# The review-assist role

## 0.1.0

The first version. The model reads the review packet of the target from the last build, acts as the critic of spec 10.4, and writes the packet with its concerns to reports/review/TAG.md with the tool write_packet. Each concern names a binding of the packet, a class of misformalization of spec 10.5 and a reason. The packet has no reviews, thus a blind re-review can use it. The run succeeds when the file has the packet of the build and each concern is valid. Review-assist never signs a review.
