package session_test

import (
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/replai/internal/session"
)

var _ = Describe("Store", func() {
	var store *session.Store

	BeforeEach(func() {
		store = session.NewStore(GinkgoT().TempDir())
	})

	It("creates sessions with unique ids", func() {
		a, err := store.Create("/proj")
		Expect(err).NotTo(HaveOccurred())
		b, err := store.Create("/proj")
		Expect(err).NotTo(HaveOccurred())
		Expect(a.ID).NotTo(Equal(b.ID))
		Expect(a.ID).To(HavePrefix("s"))
	})

	It("round-trips state through save and load", func() {
		st, err := store.Create("/proj")
		Expect(err).NotTo(HaveOccurred())
		st.Entries = append(st.Entries, &session.Entry{Kind: "stmt", Source: "x := 1", Defined: map[string]string{"x": "int"}})
		Expect(store.Save(st)).To(Succeed())

		loaded, err := store.Load(st.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(loaded.Entries).To(HaveLen(1))
		Expect(loaded.Entries[0].Source).To(Equal("x := 1"))
	})

	It("returns a helpful error for unknown sessions", func() {
		_, err := store.Load("snope")
		Expect(err).To(MatchError(ContainSubstring("replai session start")))
	})

	It("mutates state under WithLock and persists the result", func() {
		st, err := store.Create("/proj")
		Expect(err).NotTo(HaveOccurred())
		Expect(store.WithLock(st.ID, func(s *session.State) error {
			s.Entries = append(s.Entries, &session.Entry{Kind: "stmt", Source: "y := 2"})
			return nil
		})).To(Succeed())

		loaded, err := store.Load(st.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(loaded.Entries).To(HaveLen(1))
	})

	It("appends and reads log records", func() {
		st, err := store.Create("/proj")
		Expect(err).NotTo(HaveOccurred())
		rec := &session.LogRecord{Time: time.Now().UTC(), Input: "1+1", Output: json.RawMessage(`{"ok":true}`)}
		Expect(store.AppendLog(st.ID, rec)).To(Succeed())
		Expect(store.AppendLog(st.ID, rec)).To(Succeed())

		records, err := store.ReadLog(st.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(records).To(HaveLen(2))
	})

	It("deletes session files", func() {
		st, err := store.Create("/proj")
		Expect(err).NotTo(HaveOccurred())
		Expect(store.Delete(st.ID)).To(Succeed())
		_, err = store.Load(st.ID)
		Expect(err).To(HaveOccurred())
	})

	It("reports deleting an unknown session", func() {
		Expect(store.Delete("snope")).To(MatchError(ContainSubstring("not found")))
	})
})
